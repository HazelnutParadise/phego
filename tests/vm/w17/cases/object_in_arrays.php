<?php
class Counter
{
    public $n = 0;
    public $tags = [];
}

$list = [];
$list[] = new Counter;
$list[] = new Counter;
$list["named"] = new Counter;
$list[0]->n = 5;
$list[1]->n++;
$list["named"]->n += 3;
$list["named"]->tags[] = "t";
$list["named"]->tags["k"] = "v";
foreach ($list as $k => $c) {
    echo $k, ": ", $c->n, "\n";
}
echo $list["named"]->tags[0], $list["named"]->tags["k"], "\n";

$grid = [[new Counter]];
$grid[0][0]->n = 42;
$grid[0][] = new Counter;
$grid[0][1]->n = $grid[0][0]->n + 1;
echo $grid[0][0]->n, " ", $grid[0][1]->n, "\n";

$c = new Counter;
$c->tags = ["a" => new Counter];
$c->tags["a"]->n = 9;
$c->tags["a"]->tags[] = new Counter;
$c->tags["a"]->tags[0]->n = 10;
echo $c->tags["a"]->n, " ", $c->tags["a"]->tags[0]->n, "\n";

[$first, $second] = [new Counter, new Counter];
$first->n = 1;
$second->n = 2;
echo $first->n, $second->n, "\n";
['x' => $x] = ['x' => $first];
$x->n = 100;
echo "destructured handle: ", $first->n, "\n";

$same = [$first, $first];
$same[0]->n = 7;
echo "shared: ", $same[1]->n, "\n";
echo "isset element prop: ", (isset($list[0]->n) ? "yes" : "no"), (isset($list[9]->n) ? "yes" : "no"), "\n";
echo "array === with same handles: ", ([$first] === [$first] ? "yes" : "no"), "\n";
echo "array == with equal objects: ", ([new Counter] == [new Counter] ? "yes" : "no"), "\n";
echo "array === with equal objects: ", ([new Counter] === [new Counter] ? "yes" : "no"), "\n";
