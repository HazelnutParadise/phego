<?php
class P { function __call($n, $a) { echo "call $n\n"; } }
class C extends P { function __construct() { parent::__construct(); } }
new C;
