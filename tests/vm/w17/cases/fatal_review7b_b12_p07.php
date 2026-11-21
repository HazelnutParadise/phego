<?php
class P { }
class C extends P { function __construct() { $m = "__construct"; parent::$m(); } }
new C;
