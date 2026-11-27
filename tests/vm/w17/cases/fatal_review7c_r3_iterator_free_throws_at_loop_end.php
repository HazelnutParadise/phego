<?php
class T { function __destruct() { echo "T dtor\n"; throw new Exception("from iter"); } }
function f() { foreach ([new T, 2] as $v) { $v = null; try { return 1; } catch (Exception $e) { echo "caught ", $e->getMessage(), "\n"; } } echo "after loop\n"; return 9; }
echo f(), "\n";
