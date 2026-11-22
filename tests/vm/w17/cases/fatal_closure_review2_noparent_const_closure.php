<?php
class A { function t() { return function () { return parent::X; }; } }
echo "before\n";
(new A)->t()();
