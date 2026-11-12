<?php
$name = "World";
$n = 3;
echo <<<EOT
Hello $name!
Count: {$n} items
Escapes: \t tab \x41
EOT;
echo "\n";
echo <<<'RAW'
Raw $name {$n} \t
RAW;
echo "\n";
echo <<<"QUOTED"
  Quoted $name
  QUOTED;
echo "|\n";
echo <<<EOT
    indented
      more
    back
    EOT;
echo "|\n";
$empty = <<<EOT
EOT;
echo "[", $empty, "]\n";
