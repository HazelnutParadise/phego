<?php
class User
{
    public function __construct(
        public string $name,
        protected int $age = 30,
        private array $tags = [],
        public readonly string $id = "u0",
        $plain = "not promoted",
    ) {
        echo "body sees ", $this->name, " ", $plain, "\n";
        $this->name = $this->name . "!";
    }

    public function age()
    {
        return $this->age;
    }

    public function tags()
    {
        $out = "";
        foreach ($this->tags as $t) {
            $out .= "[" . $t . "]";
        }
        return $out;
    }
}

$u = new User("ann");
echo $u->name, " ", $u->age(), " '", $u->tags(), "' ", $u->id, "\n";
$u = new User("bob", "41", ["x", "y"], "u7");
echo $u->name, " ", $u->age(), " '", $u->tags(), "' ", $u->id, "\n";
$u = new User(id: "named", name: "cy");
echo $u->name, " ", $u->id, "\n";
echo "plain is not a property: ", (isset($u->plain) ? "yes" : "no"), "\n";

class Child extends User
{
    public function __construct(public $extra = "extra")
    {
        parent::__construct("child", 5);
    }
}
$c = new Child;
echo $c->name, " ", $c->age(), " ", $c->extra, " ", $c->id, "\n";

class Point
{
    public function __construct(public int $x = 0, public ?int $y = null)
    {
    }
}
$p = new Point(3);
echo $p->x, " ", ($p->y === null ? "null" : $p->y), "\n";
$p = new Point(y: 4);
echo $p->x, " ", $p->y, "\n";
foreach (new Point(1, 2) as $k => $v) {
    echo "prop ", $k, "=", $v, "\n";
}
