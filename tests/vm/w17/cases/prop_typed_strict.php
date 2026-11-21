<?php
declare(strict_types=1);

class Typed
{
    public int $i = 0;
    public float $f = 0.0;
    public ?string $s = null;
    public int|float $n = 0;
}

$t = new Typed;
$t->i = 5;
$t->f = 5;
echo "int widens to float in strict mode: ", $t->f, "\n";
$t->s = "ok";
$t->s = null;
echo "nullable null: ", ($t->s === null ? "null" : "set"), "\n";
$t->n = 1.5;
$t->n = 2;
echo "union: ", $t->n, "\n";
$t->i += 3;
echo "compound int: ", $t->i, "\n";
$t->i++;
echo "increment: ", $t->i, "\n";
$t->f *= 2;
echo "float compound: ", $t->f, "\n";
