<?php
class P
{
    public function __construct(public $a = 1, public $b = 2)
    {
    }
}
class Q
{
    public function __construct(public $a = 1, public $b = 2)
    {
    }
}
class R extends P
{
}

function show($label, $v)
{
    echo $label, ": ", ($v === true ? "true" : ($v === false ? "false" : $v)), "\n";
}

$x = new P;
$y = new P;
$z = $x;
show("same props ==", $x == $y);
show("same props ===", $x === $y);
show("same handle ===", $x === $z);
show("!= ", $x != $y);
show("!==", $x !== $y);
$y->b = 3;
show("diff props ==", $x == $y);
show("<=> less", $x <=> $y);
show("<=> greater", $y <=> $x);
show("<=> equal", $x <=> $z);
show("<", $x < $y);
show(">", $x > $y);
$q = new Q;
show("different class ==", $x == $q);
show("different class <=>", $x <=> $q);
show("subclass ==", new P == new R);
show("loose string vs numeric prop", new P("1") == new P(1));
show("loose 'abc' vs 0", new P("abc") == new P(0));
show("loose null vs false", new P(null) == new P(false));

$n1 = new P(new P(1, 2), [1, 2]);
$n2 = new P(new P(1, 2), [1, 2]);
show("nested ==", $n1 == $n2);
$n2->a->b = 5;
show("nested changed ==", $n1 == $n2);
show("nested <=>", $n1 <=> $n2);

show("object == true", $x == true);
show("object == null", $x == null);
show("object === null", $x === null);
show("object != false", $x != false);

class Dyn
{
}
$d1 = new Dyn;
$d2 = new Dyn;
show("empty objects ==", $d1 == $d2);
show("empty objects <=>", $d1 <=> $d2);
