--TEST--
WeakReference and WeakMap observe objects without owning them
--FILE--
<?php
class W { function __destruct(){ global $wr; echo "destruct sees ", ($wr->get() === null ? "null" : "object"), "\n"; } }
$o = new W();
$wr = WeakReference::create($o);
var_dump($wr === WeakReference::create($o));
var_dump($wr->get() === $o);
$map = new WeakMap(); $map[$o] = 'payload'; echo count($map), "\n";
unset($o);
var_dump($wr->get());
echo count($map), "\n";
--EXPECT--
bool(true)
bool(true)
1
destruct sees object
NULL
0
