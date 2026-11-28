<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
class C { function __construct(...$a) { echo "C ctor\n"; } function __destruct() { echo "~C\n"; } }
class K { function m(...$a) {} function __destruct() { echo "~K\n"; } }
function t() { echo "t\n"; throw new Exception("x"); }
function h1() { $c = new C(new D(1), t()); }
try { h1(); } catch (Exception $e) { echo "caught1\n"; }
function h2() { (new K)->m(new D(2), t()); }
try { h2(); } catch (Exception $e) { echo "caught2\n"; }
function h3() { $x = [new D(3), (new K)->m(new D(4), new C(new D(5), t()))]; }
try { h3(); } catch (Exception $e) { echo "caught3\n"; }
