golden.txt holds `name|flags|sha256` lines: the SVG produced by the CLI on
examples/owl.png with `-n 15 -j 1 -seed 5` plus the flags. A change here means
the output of the algorithm changed. Regenerate only when that is intended:
build the CLI, run each case, and write the new sha256 of the SVG.
