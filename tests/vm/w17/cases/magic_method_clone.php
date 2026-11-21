<?php
class Address
{
    public function __construct(public $city)
    {
    }
}

class Person
{
    public $tags = ["a"];
    public static $clones = 0;

    public function __construct(public $name, public Address $addr)
    {
    }
}

class DeepPerson extends Person
{
    public function __clone()
    {
        echo "__clone on ", $this->name, "\n";
        $this->addr = clone $this->addr;
        $this->name .= " (copy)";
        static::$clones++;
    }
}

$p = new Person("ann", new Address("Paris"));
$q = clone $p;
$q->name = "bob";
$q->tags[] = "b";
$q->addr->city = "Rome";
echo "shallow: ", $p->name, " ", $p->addr->city, " tags ", $p->tags[0], (isset($p->tags[1]) ? "+" : ""), "\n";
echo "same address object: ", ($p->addr === $q->addr ? "yes" : "no"), "\n";
echo "equal but not identical: ", ($p == clone $p ? "==" : "!="), " ", ($p === clone $p ? "===" : "!=="), "\n";

$d = new DeepPerson("cy", new Address("Oslo"));
$e = clone $d;
$e->addr->city = "Bergen";
echo "deep: ", $d->name, " ", $d->addr->city, " | ", $e->name, " ", $e->addr->city, "\n";
$f = clone $e;
echo "clone count: ", DeepPerson::$clones, " ", $f->name, "\n";

$p->dyn = "dynamic";
$g = clone $p;
echo "dynamic props copied: ", $g->dyn, "\n";
echo "clone of new: ", (clone new Address("Lima"))->city, "\n";
echo "class kept: ", (clone $d)::class, "\n";
