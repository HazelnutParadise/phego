<?php
class A implements IteratorAggregate { function getIterator() { return [1, 2]; } }
try { foreach (new A as $v) {} } catch (Exception $e) { echo $e::class, ": ", $e->getMessage(), "\n"; }
