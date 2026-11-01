--TEST--
Destructor runs after the last object reference is removed
--FILE--
<?php
class W06Object {
    public function __destruct() { echo "destruct\n"; }
}
$first = new W06Object();
$second = $first;
unset($first);
echo "after first\n";
unset($second);
echo "after second\n";
?>
--EXPECT--
after first
destruct
after second
