--TEST--
Known environment skip
--SKIPIF--
<?php echo "skip synthetic unavailable environment"; ?>
--FILE--
<?php echo "unreachable\n"; ?>
--EXPECT--
unreachable
