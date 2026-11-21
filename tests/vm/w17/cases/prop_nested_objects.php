<?php
class Node
{
    public $child;
    public $value = 0;
    public $items = [];
}

$root = new Node;
$root->child = new Node;
$root->child->child = new Node;
$root->child->child->value = 3;
$root->child->value = 2;
$root->value = 1;
echo $root->value, $root->child->value, $root->child->child->value, "\n";
$root->child->child->value += 10;
$root->child->value++;
--$root->value;
echo $root->value, " ", $root->child->value, " ", $root->child->child->value, "\n";
$root->child->items[] = "a";
$root->child->items["x"]["y"] = "b";
echo $root->child->items[0], $root->child->items["x"]["y"], "\n";
$root->child->items[0] .= "!";
echo $root->child->items[0], "\n";

$alias = $root->child;
$alias->value = 99;
echo "alias: ", $root->child->value, "\n";
$root->child = null;
echo "alias survives: ", $alias->value, " ", $alias->child->value, "\n";

$root->items["obj"] = new Node;
$root->items["obj"]->value = "in array in object";
echo $root->items["obj"]->value, "\n";
echo "isset deep: ", (isset($alias->child->value) ? "yes" : "no"), (isset($root->child->value) ? "yes" : "no"), "\n";
echo "coalesce deep: ", ($root->child->child->value ?? "none"), "\n";
