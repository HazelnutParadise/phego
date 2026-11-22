<?php
function f() {
  throw
    new
      LogicException("l");
}
try { f(); } catch (Exception $e) { echo $e->getLine(), "\n"; }
