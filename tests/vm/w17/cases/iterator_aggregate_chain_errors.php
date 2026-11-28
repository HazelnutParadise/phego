<?php
class A implements IteratorAggregate { #[\ReturnTypeWillChange] function getIterator() { return new B; } }
class B implements IteratorAggregate { #[\ReturnTypeWillChange] function getIterator() { return new stdClass; } }
try { foreach (new A as $v) {} } catch (Exception $e) { echo $e::class, ": ", $e->getMessage(), "\n"; }
class C implements IteratorAggregate { function getIterator(): Traversable { throw new LogicException("no"); } }
try { foreach (new C as $v) {} } catch (LogicException $e) { echo "caught ", $e->getMessage(), "\n"; }
