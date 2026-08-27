# Brand assets

`icon-source.jpeg` is the generated sheet the icons came from: an app tile, plus
sample favicons and alternate layouts.

`app-icon-512.png` is that tile, cropped and downscaled — the icon to use where
there is room for it (README, a GitHub social preview, a PWA manifest).

`../../ui/public/favicon.svg` is **not** a downscale of it, deliberately. At 16px
the illustration's monitor, pod network, palm and sandcastle collapse into a
teal-and-tan smudge, so the favicon is a simplified mark that keeps the three
things which still read at that size: the teal tile, the sand, and one pod. The
colours are sampled from the illustration.

`../../ui/public/favicon.ico` holds 16/32/48 rasters of that mark for browsers
that will not take an SVG. Its rounded corners are filled with the frame teal
rather than left transparent, because an ICO of this form has no alpha and white
corners show as notches on a dark tab strip.
