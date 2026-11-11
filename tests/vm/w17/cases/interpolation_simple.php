<?php
$x = "val";
$i = 7;
$f = 1.5;
$b = true;
$z = null;
echo "plain $x end\n";
echo "braced {$x}end\n";
echo "int $i float $f bool $b null [$z]\n";
echo "adjacent $x$i\n";
echo "not a var: $ 5 and \$x\n";
echo "brace text { $x }\n";
