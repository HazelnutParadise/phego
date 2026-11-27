<?php
$a = null;
try {
  throw $a
    ??
    5;
} catch (Error $e) { echo $e->getMessage(), " ", $e->getLine(), "\n"; }
