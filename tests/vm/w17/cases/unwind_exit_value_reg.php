<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
$x = [new D(1), exit("bye\n")];
