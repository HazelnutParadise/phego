<?php
abstract class X implements Iterator {}
class Y extends X implements IteratorAggregate { function getIterator(): Iterator { return $this; } }
