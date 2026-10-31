--TEST--
A destructor exception from the iterated array's final release escapes foreach
--FILE--
<?php
class T { function __destruct() { throw new Exception("from destructor"); } }
$list = [new T, 1];
try { foreach ($list as &$item) { unset($list); } echo "no exception\n"; } catch (Exception $e) { echo "caught ", $e->getMessage(), "\n"; }
--EXPECT--
caught from destructor
