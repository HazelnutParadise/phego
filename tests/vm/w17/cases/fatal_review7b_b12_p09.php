<?php
class A { function f() { static::__construct(); } }
(new A)->f();
