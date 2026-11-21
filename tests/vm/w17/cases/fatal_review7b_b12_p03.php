<?php
class A { function f() { A::__construct(); } }
(new A)->f();
