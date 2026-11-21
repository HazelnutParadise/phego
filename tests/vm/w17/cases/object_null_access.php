<?php
$n = null;
echo "read prop on null: [", $n->prop, "]\n";
$i = 5;
echo "read prop on int: [", $i->prop, "]\n";
$s = "str";
echo "read prop on string: [", $s->prop, "]\n";
$a = [1];
echo "read prop on array: [", $a->prop, "]\n";
$f = false;
echo "read prop on false: [", $f->prop, "]\n";
echo "read prop on undefined: [", $undef->prop, "]\n";
echo "isset on null: ", (isset($n->prop) ? "yes" : "no"), "\n";
echo "isset nested: ", (isset($n->a->b->c) ? "yes" : "no"), "\n";
echo "empty on null: ", (empty($n->prop) ? "yes" : "no"), "\n";
echo "coalesce: ", ($n->prop ?? "default"), "\n";
echo "coalesce nested: ", ($n->a->b ?? "deep default"), "\n";
unset($n->prop);
echo "unset on null is silent\n";

class Holder
{
    public $inner = null;
}
$h = new Holder;
echo "chain through null property: [", $h->inner->x, "]\n";
echo "isset chain: ", (isset($h->inner->x) ? "yes" : "no"), "\n";
echo "nullsafe: [", $h->inner?->x, "]\n";
function f()
{
    $local = null;
    return $local->field;
}
echo "in function: [", f(), "]\n";
