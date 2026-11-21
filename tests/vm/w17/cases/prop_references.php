<?php
class Holder
{
    public $v = 1;
    public $arr = [1, 2, 3];
    public int $typed = 5;
}

function add_one(&$x)
{
    $x++;
}

function append(&$list, $v)
{
    $list[] = $v;
}

$h = new Holder;
$r = &$h->v;
$r = 10;
echo "write through reference: ", $h->v, "\n";
$h->v = 20;
echo "read through reference: ", $r, "\n";
unset($r);
$h->v = 30;
echo "after unset ref: ", $h->v, "\n";

add_one($h->v);
echo "by-ref param: ", $h->v, "\n";
append($h->arr, 4);
echo "by-ref array param: ", $h->arr[3], "\n";
add_one($h->arr[0]);
echo "by-ref element: ", $h->arr[0], "\n";

foreach ($h->arr as &$item) {
    $item *= 10;
}
unset($item);
echo "foreach by ref: ", $h->arr[0], " ", $h->arr[3], "\n";

$t = &$h->typed;
$t = "8";
echo "typed reference coerces: ", $h->typed, "\n";
add_one($h->typed);
echo "typed by-ref param: ", $h->typed, "\n";

$a = new Holder;
$b = new Holder;
$a->v = &$b->v;
$b->v = "shared";
echo "property to property ref: ", $a->v, "\n";
$copy = clone $a;
$copy->v = "copy writes the shared reference";
echo "clone keeps reference: ", $b->v, "\n";

$local = "local";
$h->v = &$local;
$local = "changed local";
echo "property bound to local: ", $h->v, "\n";

$created = new Holder;
$ref = &$created->newDyn;
$ref = "via ref";
echo "reference creates dynamic: ", $created->newDyn, "\n";
add_one($created->counter);
echo "by-ref creates: ", $created->counter, "\n";
