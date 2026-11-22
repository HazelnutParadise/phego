<?php
$e = null;
try {
  throw
    $e;
} catch (Error $x) { echo $x->getLine(), "\n"; }
