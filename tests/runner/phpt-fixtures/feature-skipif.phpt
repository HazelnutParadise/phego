--TEST--
Feature precondition for classification
--SKIPIF--
<?php if (!function_exists('strlen')) echo 'skip strlen unavailable'; ?>
--FILE--
<?php echo strlen('abc'), "\n"; ?>
--EXPECT--
3
