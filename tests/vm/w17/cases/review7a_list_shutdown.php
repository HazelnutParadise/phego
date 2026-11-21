<?php
class N { public $next; function __construct(public $n) {} function __destruct() { echo "~{$this->n}\n"; } }
$head = new N(1);
$head->next = new N(2);
$tail = $head->next;
echo "end\n";
