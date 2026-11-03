--TEST--
Unexpected pass for expected failure
--XFAIL--
Synthetic expected mismatch
--FILE--
<?php echo "matching\n"; ?>
--EXPECT--
matching
