<?php
interface I { const X = 1; }
class A { private const X = 2; }
class B extends A implements I { }
echo B::X, "\n";
