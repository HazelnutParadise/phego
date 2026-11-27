<?php
function mk($m) { return $m; }
try {
  throw mk(
    "s"
  );
} catch (Error $e) { echo $e->getMessage(), " ", $e->getLine(), "\n"; }
