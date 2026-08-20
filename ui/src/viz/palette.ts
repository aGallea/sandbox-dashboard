/**
 * The dashboard's chart tokens, in one place so every page draws from the same
 * system.
 *
 * Validated with the data-viz palette validator against the #ffffff card
 * surface: the five categorical slots pass every gate (worst adjacent CVD
 * ΔE 9.1, normal-vision ΔE 19.6), as does the five-step ordinal blue ramp.
 * Three of the categorical fills sit under 3:1 on white, so a chart using them
 * ships visible labels — a legend with values, or labels on the marks.
 *
 * Assign the categorical slots in this order and never cycle past the last one:
 * a sixth identity folds into Other instead of inventing a hue.
 */
export const SERIES = ['#2a78d6', '#eb6834', '#1baf7a', '#eda100', '#e87ba4'];

/** The folded tail is a leftover, not an identity — it never takes a hue. */
export const OTHER_COLOR = '#94a3b8';

/** For ordered categories: one hue, light to dark, rather than five identities. */
export const AGE_RAMP = ['#86b6ef', '#5598e7', '#2a78d6', '#1c5cab', '#104281'];

/**
 * Reserved for state, never for a series. Each is paired with an icon and a
 * word wherever it appears, so meaning never rests on hue alone.
 */
export const STATUS = {
  ready: '#0ca30c',
  pending: '#fab219',
  failed: '#d03b3b',
  idle: '#94a3b8',
};

/** Chart chrome: recessive hairlines, one shade off the surface. */
export const AXIS = '#94a3b8';
export const GRID = '#e2e8f0';
export const HOVER = '#f8fafc';

export const tooltipStyle = {
  fontSize: 12,
  borderRadius: 8,
  border: `1px solid ${GRID}`,
  boxShadow: 'none',
  padding: '4px 8px',
};
