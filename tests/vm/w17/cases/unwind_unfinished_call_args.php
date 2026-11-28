<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
class K { function m(...$a) { echo "m\n"; } }
class C { function __construct(...$a) {} }
function t() { echo "t\n"; throw new Exception("x"); }
function g(...$a) { return 1; }
function h1() { (new K)->m(new D(1), t()); }
try { h1(); } catch (Exception $e) { echo "caught1\n"; }
function h2() { new C(new D(2), t()); }
try { h2(); } catch (Exception $e) { echo "caught2\n"; }
function h3() { g(new D(3), g(new D(4), t())); }
try { h3(); } catch (Exception $e) { echo "caught3\n"; }
try { $x = [new D(5), g(new D(6), t())]; } catch (Exception $e) { echo "caught4\n"; }
function h5() { g(a: new D(7), b: t()); }
try { h5(); } catch (Exception $e) { echo "caught5\n"; }
function h6() { (new K)->m(new D(8), t()); }
function h6o() { try { h6(); } finally { echo "fin6\n"; } }
try { h6o(); } catch (Exception $e) { echo "caught6 ", $e->getTraceAsString(), "\n"; }
