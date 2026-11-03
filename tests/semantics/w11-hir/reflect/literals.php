<?php
$values[] = 0x7FFF_FFFF;
$values[] = 0b1010;
$values[] = 0o17;
$values[] = 017;
$values[] = 1_000_000;
$values[] = 1.5e3;
$values[] = .5;
$values[] = 1e400;
$values[] = 0x1FFFFFFFFFFFFFFFF;
$values[] = 0b11111111111111111111111111111111111111111111111111111111111111111;
$values[] = 0777777777777777777777777;
$values[] = 9223372036854775807;
$values[] = 9223372036854775808;
$values[] = 'single \' \\ \n $x';
$values[] = "double \t\n\r\v\e\f\\\$\"";
$values[] = "hex \x41\x4 \xZ oct \101\7\400 uni \u{1F600}\u{48} \u not";
$values[] = "keep \q \' \`";
$values[] = <<<EOT
    indented
      more
    last
    EOT;
$values[] = <<<"EOT"
  a\tb
  "quoted" \"
  EOT;
$values[] = <<<'EOT'
    raw \t $x
     kept
    EOT;
$values[] = <<<EOT
EOT;
$values[] = <<<EOT
  x

  y
  EOT;
$values[] = b"bin\n";
$values[] = true;
$values[] = NULL;
$values[] = False;
