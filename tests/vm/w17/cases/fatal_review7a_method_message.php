<?php
class A { function m(int $a, string ...$rest) {} }
(new A)->m(1, "a", []);
