<?php
class Res
{
    public function __construct(public $name)
    {
        echo "open ", $name, "\n";
    }

    public function __destruct()
    {
        echo "close ", $this->name, "\n";
    }
}

$a = new Res("a");
unset($a);
echo "after unset a\n";

$b = new Res("b");
$b = new Res("b2");
echo "after reassign b\n";

$c = new Res("c");
$c = null;
echo "after null c\n";

$d = new Res("d");
$d2 = $d;
unset($d);
echo "d still referenced\n";
$d2 = 1;
echo "after last handle of d\n";

$e = new Res("e");
$ref = &$e;
unset($e);
echo "e alive through reference\n";
unset($ref);
echo "after reference gone\n";

$list = [new Res("x"), new Res("y")];
unset($list[0]);
echo "after unset element\n";
$list = [];
echo "after array cleared\n";

$o = new Res("holder");
$o->child = null;
echo "end of script\n";
