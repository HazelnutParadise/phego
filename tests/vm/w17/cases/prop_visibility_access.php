<?php
class Account
{
    public $owner = "ann";
    protected $balance = 100;
    private $pin = "1234";

    public function summary()
    {
        return $this->owner . " " . $this->balance . " " . $this->pin;
    }

    public function peekOther(Account $o)
    {
        return "other pin " . $o->pin . " balance " . $o->balance;
    }

    public function setPinOn(Account $o, $pin)
    {
        $o->pin = $pin;
    }
}

class Savings extends Account
{
    public function addInterest()
    {
        $this->balance += 5;
        return $this->balance;
    }

    public function siblingBalance(Checking $c)
    {
        return "sibling balance " . $c->balance;
    }
}

class Checking extends Account
{
}

$a = new Account;
$b = new Account;
echo $a->summary(), "\n";
echo $a->peekOther($b), "\n";
$a->setPinOn($b, "9999");
echo $b->summary(), "\n";
$s = new Savings;
echo "subclass protected: ", $s->addInterest(), "\n";
echo "sibling via common ancestor: ", $s->siblingBalance(new Checking), "\n";
echo "base method on subclass: ", $s->summary(), "\n";
echo "isset from outside: ", (isset($a->balance) ? "yes" : "no"), (isset($a->pin) ? "yes" : "no"), (isset($a->owner) ? "yes" : "no"), "\n";
echo "empty from outside: ", (empty($a->balance) ? "yes" : "no"), "\n";
echo "coalesce from outside: ", ($a->pin ?? "hidden"), "\n";
