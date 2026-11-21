<?php
class Base
{
    public $pub = "p";
    protected $prot = "q";
    private $priv = "r";
    public $nested = [1, 2];
}
class Child extends Base
{
    private $priv = "child r";
    public $extra = null;
}

$o = new Child;
$o->dyn = "dynamic";
$a = (array)$o;
$n = 0;
foreach ($a as $k => $v) {
    $n++;
    echo "[", $k, "] => ", (is_array_like($v) ? "array" : $v), "\n";
}
echo "entries: ", $n, "\n";
echo "public key readable: ", $a["pub"], " ", $a["dyn"], "\n";
echo "mangled protected key: ", $a["\0*\0prot"], "\n";
echo "mangled private keys: ", $a["\0Base\0priv"], " / ", $a["\0Child\0priv"], "\n";
echo "plain private key missing: ", (isset($a["priv"]) ? "yes" : "no"), "\n";

$a["pub"] = "changed in array";
echo "array is a copy: ", $o->pub, "\n";
$o->nested[] = 3;
echo "copy keeps old nested: ", $a["nested"][1], " ", (isset($a["nested"][2]) ? "has 3" : "no 3"), "\n";

function is_array_like($v)
{
    return $v === [1, 2];
}

$empty = (array)new class_with_nothing;
echo "empty cast is empty: ", ($empty === [] ? "yes" : "no"), "\n";
class class_with_nothing
{
}

$arr = [1, 2];
echo "array cast of array is identity: ", ((array)$arr === $arr ? "yes" : "no"), "\n";
$s = (array)"str";
echo "scalar cast: ", $s[0], "\n";
$nul = (array)null;
echo "null cast is empty: ", ($nul === [] ? "yes" : "no"), "\n";
