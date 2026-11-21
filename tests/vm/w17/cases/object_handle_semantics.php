<?php
class Item
{
    public $v = 1;
    public $list = [];
}

function mutate($o)
{
    $o->v = 100;
    $o->list[] = "added";
}

function reassign($o)
{
    $o = new Item;
    $o->v = -1;
    return $o;
}

function reassign_by_ref(&$o)
{
    $o = new Item;
    $o->v = 55;
}

$a = new Item;
mutate($a);
echo "after mutate: ", $a->v, " ", $a->list[0], "\n";
$r = reassign($a);
echo "after reassign: ", $a->v, " returned: ", $r->v, "\n";
reassign_by_ref($a);
echo "after by-ref reassign: ", $a->v, "\n";

$b = $a;
$b->v = 7;
echo "alias: ", $a->v, " same: ", ($a === $b ? "yes" : "no"), "\n";
$b = new Item;
echo "rebinding b leaves a: ", $a->v, " ", $b->v, "\n";

$c = &$a;
$c = new Item;
echo "reference rebinding changes a: ", $a->v, "\n";

$arr = [$a, $a];
$arr[0]->v = 9;
echo "array holds handles: ", $arr[1]->v, " ", $a->v, "\n";
$copy = $arr;
$copy[0]->v = 10;
echo "array copy still shares objects: ", $arr[0]->v, "\n";

$list = [new Item, new Item];
foreach ($list as $item) {
    $item->v++;
}
echo "foreach by value mutates objects: ", $list[0]->v, $list[1]->v, "\n";
foreach ($list as $item) {
    $item = new Item;
    $item->v = 99;
}
echo "foreach rebinding does not: ", $list[0]->v, "\n";

class Node
{
    public $next = null;
    public function __construct(public $name)
    {
    }
}
$first = new Node("one");
$first->next = new Node("two");
$first->next->next = new Node("three");
$n = $first;
$names = "";
while ($n !== null) {
    $names .= $n->name . ">";
    $n = $n->next;
}
echo $names, "\n";
$first->next->next->next = $first;
echo "cycle: ", $first->next->next->next->next->name, "\n";
