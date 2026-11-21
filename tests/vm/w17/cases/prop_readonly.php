<?php
class Money
{
    public function __construct(
        public readonly int $amount,
        public readonly string $currency = "EUR",
    ) {
    }

    public function withAmount($a)
    {
        return new static($a, $this->currency);
    }
}

class Lazy
{
    public readonly array $items;
    public readonly ?string $late;

    public function init()
    {
        $this->items = [1, 2];
        return $this;
    }

    public function isReady()
    {
        return isset($this->items) ? "ready" : "not ready";
    }

    public function setLate($v)
    {
        $this->late = $v;
    }
}

class Cloned
{
    public function __construct(public readonly int $v, public readonly array $list = [])
    {
    }

    public function __clone()
    {
        $this->v = $this->v + 100;
    }
}

$m = new Money(5);
echo $m->amount, " ", $m->currency, "\n";
$m2 = $m->withAmount(10);
echo $m2->amount, " ", $m2->currency, " original ", $m->amount, "\n";

$l = new Lazy;
echo $l->isReady(), "\n";
echo $l->init()->isReady(), " ", $l->items[1], "\n";
echo "uninitialized readonly isset: ", (isset($l->late) ? "yes" : "no"), "\n";
$l->setLate(null);
echo "initialized to null isset: ", (isset($l->late) ? "yes" : "no"), "\n";

$c = new Cloned(1, ["a"]);
$d = clone $c;
echo "clone reinitialized in __clone: ", $c->v, " ", $d->v, " ", $d->list[0], "\n";

$obj = new Cloned(5, [new Money(1)]);
echo "inner object of readonly array is mutable via handle: ", $obj->list[0]->amount, "\n";
$copy = $obj->list;
$copy[] = "x";
echo "copy of readonly array is free: ", (isset($copy[1]) ? "yes" : "no"), "\n";
foreach ($m as $k => $v) {
    echo "iterate ", $k, "=", $v, "\n";
}
