<?php
class C { function __destruct() { echo "~C\n"; } }
class D { public $c; function __construct() { $this->c = new C; $u = null; $u->x(); } function __destruct() { echo "~D\n"; } }
$d = new D;
