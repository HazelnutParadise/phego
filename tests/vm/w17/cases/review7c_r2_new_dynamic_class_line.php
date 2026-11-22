<?php
$x = null;
$e = new ($x
  ?? "Exception")("m");
echo $e->getLine(), "\n";
