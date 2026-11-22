<?php
// CVE: CVE-2010-4697
// The object on which __get, __set, __isset or __unset ran could be freed
// while the handler still used it, when the only reference was overwritten
// inside the handler (Zend/tests/magic_methods/bug52879.phpt).
class MyClass {
    public $myRef;
    public function __set($property, $value) {
        $this->myRef = $value;
    }
}
$myGlobal = new MyClass();
$myGlobal->myRef = &$myGlobal;
$myGlobal->myNonExistentProperty = "ok\n";
echo $myGlobal;

class Getter {
    public $self;
    public function __get($name) {
        $this->self = "replaced";
        return $name . " read";
    }
    public function __isset($name) {
        $this->self = null;
        return true;
    }
    public function __unset($name) {
        $this->self = 0;
        echo "unset ", $name, "\n";
    }
}
$g = new Getter();
$g->self = &$g;
echo $g->missing, "\n";
echo $g, "\n";
$h = new Getter();
$h->self = &$h;
echo isset($h->missing) ? "isset\n" : "not set\n";
var_dump_null($h);
$u = new Getter();
$u->self = &$u;
unset($u->missing);
echo $u, "\n";
function var_dump_null($x) {
    echo $x === null ? "null\n" : "not null\n";
}
