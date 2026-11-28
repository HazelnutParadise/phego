<?php
class A implements IteratorAggregate { function getIterator(): Iterator { return (function () { yield 1; })(); } }
foreach (new A as &$v) {}
