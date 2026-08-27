import { describe, expect, it } from 'vitest';

import { guessLevel, normaliseLevel, parseLine, parseLogs, shortTime } from './format';

describe('parseLine', () => {
  it('renders a slog-style JSON line as time, level, message and the rest as k=v', () => {
    const line = parseLine(
      '{"time":"2026-08-27T10:11:12.345Z","level":"ERROR","msg":"pull failed","image":"ghcr.io/x:1","attempt":3}',
    );
    expect(line.level).toBe('error');
    expect(line.text).toBe(
      `${shortTime('2026-08-27T10:11:12.345Z')} ERROR pull failed image=ghcr.io/x:1 attempt=3`,
    );
  });

  it('understands zap epoch seconds and pino numeric levels', () => {
    const zap = parseLine('{"ts":1724745600.5,"level":"warn","msg":"slow"}');
    expect(zap.level).toBe('warn');
    expect(zap.text.startsWith(shortTime(1724745600.5))).toBe(true);

    const pino = parseLine('{"level":50,"time":1724745600500,"msg":"boom"}');
    expect(pino.level).toBe('error');
    expect(pino.text).toContain('ERROR boom');
  });

  it('quotes values that would not survive as a bare token', () => {
    const line = parseLine('{"msg":"hi","path":"a b","n":{"x":1}}');
    expect(line.text).toBe('hi path="a b" n={"x":1}');
  });

  it('leaves a plain line alone and guesses its level from the text', () => {
    expect(parseLine('E0827 10:11:12.000 1 main.go:1] klog error').level).toBe('error');
    expect(parseLine('[WARN] disk almost full')).toEqual({
      raw: '[WARN] disk almost full',
      level: 'warn',
      text: '[WARN] disk almost full',
    });
    expect(parseLine('level=info msg=started').level).toBe('info');
    expect(parseLine('just some output').level).toBe('unknown');
  });

  it('keeps a line that only looks like JSON', () => {
    const raw = '{not json';
    expect(parseLine(raw)).toEqual({ raw, level: 'unknown', text: raw });
    expect(parseLine('{"a":1}').text).toBe('a=1');
  });
});

describe('normaliseLevel and guessLevel', () => {
  it('maps the common spellings onto four buckets', () => {
    expect(normaliseLevel('CRITICAL')).toBe('error');
    expect(normaliseLevel('Warning')).toBe('warn');
    expect(normaliseLevel('trace')).toBe('debug');
    expect(normaliseLevel('NOTICE')).toBe('info');
    expect(normaliseLevel('DEFAULT')).toBe('unknown');
    expect(guessLevel('W0827 x')).toBe('warn');
  });
});

describe('parseLogs', () => {
  it('drops only the trailing newline', () => {
    expect(parseLogs('a\n\nb\n').map((l) => l.raw)).toEqual(['a', '', 'b']);
    expect(parseLogs('')).toEqual([]);
  });
});
