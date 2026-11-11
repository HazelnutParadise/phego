--TEST--
Formatted output placeholder
--FILE--
<?php echo "items: 42\n"; ?>
--EXPECTF--
items: %d
