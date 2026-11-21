<?php
class C
{
    public $n = 5;
    public $s = "a";
    public $null = null;
    public $f = 1.5;
    public $arr = [1];
}

$c = new C;
echo "post inc returns old: ", $c->n++, " now ", $c->n, "\n";
echo "pre inc returns new: ", ++$c->n, "\n";
echo "post dec: ", $c->n--, " pre dec: ", --$c->n, "\n";
$c->n += 10;
$c->n -= 3;
$c->n *= 2;
$c->n /= 4;
echo "arith: ", $c->n, "\n";
$c->n = 17;
$c->n %= 5;
$c->n **= 3;
echo "mod pow: ", $c->n, "\n";
$c->n = 6;
$c->n |= 1;
$c->n &= 5;
$c->n ^= 3;
$c->n <<= 2;
$c->n >>= 1;
echo "bitwise: ", $c->n, "\n";
$c->s .= "b";
$c->s .= 3;
echo "concat: ", $c->s, "\n";
$c->s++;
echo "string increment: ", $c->s, "\n";
$c->null++;
echo "null increment: ", $c->null, "\n";
$x = new C;
$x->null--;
echo "null decrement stays null: ", ($x->null === null ? "null" : $x->null), "\n";
$c->f++;
echo "float inc: ", $c->f, "\n";
$c->null ??= "coalesced";
echo "??= on non-null: ", $c->null, "\n";
$x->null ??= "coalesced";
echo "??= on null: ", $x->null, "\n";
$x->undefinedDyn ??= "created";
echo "??= on undefined creates: ", $x->undefinedDyn, "\n";
$c->undef += 1;
echo "compound on undefined: ", $c->undef, "\n";
$c->arr += [5, 6, 7];
echo "array union: ", $c->arr[0], $c->arr[1], $c->arr[2], "\n";

function side($v)
{
    echo "[side ", $v, "]";
    return $v;
}
$c->n = 1;
$c->n += side(2);
echo " ", $c->n, "\n";
$y = new C;
$y->null ??= side("only once");
$y->null ??= side("never");
echo " ", $y->null, "\n";
