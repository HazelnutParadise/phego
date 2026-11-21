<?php
class MathX
{
    public function fact($n)
    {
        return $n <= 1 ? 1 : $n * $this->fact($n - 1);
    }

    public static function fib($n)
    {
        return $n < 2 ? $n : static::fib($n - 1) + self::fib($n - 2);
    }

    public function isEven($n)
    {
        return $n === 0 ? true : $this->isOdd($n - 1);
    }

    public function isOdd($n)
    {
        return $n === 0 ? false : $this->isEven($n - 1);
    }
}

class Tree
{
    public $left;
    public $right;
    public function __construct(public $v, $l = null, $r = null)
    {
        $this->left = $l;
        $this->right = $r;
    }
    public function sum()
    {
        return $this->v + ($this->left?->sum() ?? 0) + ($this->right?->sum() ?? 0);
    }
    public function depth()
    {
        $l = $this->left ? $this->left->depth() : 0;
        $r = $this->right ? $this->right->depth() : 0;
        return 1 + ($l > $r ? $l : $r);
    }
}

$m = new MathX;
echo $m->fact(10), "\n";
echo MathX::fib(15), "\n";
echo ($m->isEven(10) ? "even" : "odd"), " ", ($m->isOdd(7) ? "odd" : "even"), "\n";
$t = new Tree(1, new Tree(2, new Tree(4)), new Tree(3, null, new Tree(5, new Tree(6))));
echo $t->sum(), " ", $t->depth(), "\n";
