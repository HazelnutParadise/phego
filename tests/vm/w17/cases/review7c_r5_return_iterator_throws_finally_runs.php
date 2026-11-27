<?php
class T { function __destruct(){ echo "~T\n"; throw new Exception("t"); } }
function f() { foreach ([1] as $v) { try { foreach ([new T] as $w) { $w = null; return "r"; } } finally { echo "F\n"; } } }
try { echo f(), "\n"; } catch (Exception $e) { echo "G ", $e->getMessage(), "\n"; }
