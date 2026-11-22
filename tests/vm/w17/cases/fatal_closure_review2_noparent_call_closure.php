<?php
class A { function t() { return fn() => parent::m(); } }
echo "before\n";
(new A)->t()();
