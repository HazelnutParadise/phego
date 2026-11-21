<?php
class Strict
{
    public $declared = 1;
}

$s = new Strict;
$s->declared = 2;
echo "declared write is silent\n";
$s->extra = "first";
echo "dynamic value: ", $s->extra, "\n";
$s->extra = "second";
echo "second write to same dynamic property is silent: ", $s->extra, "\n";
$t = new Strict;
$t->extra = "other instance";
echo "each instance warns\n";
unset($s->declared);
$s->declared = 3;
echo "re-adding unset declared property is silent: ", $s->declared, "\n";

class Inner
{
    public function addSelf()
    {
        $this->fromInside = "inside";
        return $this->fromInside;
    }
}
echo (new Inner)->addSelf(), "\n";

function in_function($o)
{
    $o->viaFunction = 1;
}
in_function(new Strict);

$s->arr[] = "dynamic array append";
echo $s->arr[0], "\n";
$s->counter ??= 10;
echo "coalesce assign: ", $s->counter, "\n";
$r = &$s->byRef;
echo "reference creates: ", ($s->byRef === null ? "null" : "set"), "\n";
$m = "nameVar";
$s->$m = "variable name";
echo $s->nameVar, "\n";
