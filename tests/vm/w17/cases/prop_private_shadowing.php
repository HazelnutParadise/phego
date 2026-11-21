<?php
class ParentC
{
    private $secret = "parent secret";
    public $shared = "parent shared";

    public function parentSees()
    {
        return $this->secret;
    }

    public function setParentSecret($v)
    {
        $this->secret = $v;
    }
}

class ChildC extends ParentC
{
    public $secret = "child public";

    public function childSees()
    {
        return $this->secret;
    }
}

class GrandC extends ParentC
{
    public function grandReads()
    {
        return $this->secret ?? "not visible";
    }

    public function grandWrites()
    {
        $this->secret = "grand dynamic";
        return $this->secret . " / " . $this->parentSees();
    }
}

$c = new ChildC;
echo "parent method: ", $c->parentSees(), "\n";
echo "child method: ", $c->childSees(), "\n";
echo "outside: ", $c->secret, "\n";
$c->setParentSecret("changed parent copy");
echo "after parent write: ", $c->parentSees(), " | ", $c->secret, "\n";
$c->secret = "changed child copy";
echo "after outside write: ", $c->parentSees(), " | ", $c->secret, "\n";
$n = 0;
foreach ((array)$c as $k => $v) {
    $n++;
}
echo "two slots for secret plus shared: ", $n, "\n";

$g = new GrandC;
echo "grand read: ", $g->grandReads(), "\n";
echo "grand write: ", $g->grandWrites(), "\n";
