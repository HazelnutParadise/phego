<?php
class Animal
{
    protected $sound = "...";
    public $name;

    public function __construct($name)
    {
        $this->name = $name;
        echo "Animal ctor ", $name, "\n";
    }

    public function speak()
    {
        return $this->name . " says " . $this->sound;
    }

    public function kind()
    {
        return "animal";
    }
}

class Dog extends Animal
{
    protected $sound = "woof";

    public function kind()
    {
        return "dog, a kind of " . parent::kind();
    }
}

class Puppy extends Dog
{
    public function __construct($name)
    {
        echo "Puppy ctor before parent\n";
        parent::__construct("little " . $name);
    }

    public function speak()
    {
        return parent::speak() . " (squeaky)";
    }

    public function kind()
    {
        return "puppy, " . parent::kind();
    }
}

class Silent extends Animal
{
    public function __construct()
    {
        echo "Silent ctor without parent call\n";
    }
}

$d = new Dog("rex");
echo $d->speak(), "\n";
echo $d->kind(), "\n";
$p = new Puppy("bo");
echo $p->speak(), "\n";
echo $p->kind(), "\n";
$s = new Silent;
echo "name is null: ", ($s->name === null ? "yes" : "no"), "\n";
echo $s->speak(), "\n";

echo "instanceof chain: ",
    ($p instanceof Puppy ? "P" : "-"),
    ($p instanceof Dog ? "D" : "-"),
    ($p instanceof Animal ? "A" : "-"),
    ($d instanceof Puppy ? "P" : "-"), "\n";
