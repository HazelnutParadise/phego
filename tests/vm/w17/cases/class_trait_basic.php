<?php
trait Greets
{
    public $greeting = "hello";

    public function greet($who)
    {
        return $this->greeting . " " . $who . " from " . self::class . "/" . static::class;
    }
}

trait Counts
{
    private $count = 0;

    public function bump()
    {
        $this->count++;
        return $this;
    }

    public function count()
    {
        return $this->count;
    }
}

class Person
{
    use Greets, Counts;
}

class Robot
{
    use Greets;
    public $greeting = "hello";
}

class LoudPerson extends Person
{
    public function greet($who)
    {
        return "LOUD " . parent::greet($who);
    }
}

$p = new Person;
echo $p->greet("world"), "\n";
echo $p->bump()->bump()->count(), "\n";
$r = new Robot;
echo $r->greet("human"), "\n";
$l = new LoudPerson;
echo $l->greet("all"), "\n";
echo "trait method __CLASS__-like: ", $l->bump()->count(), "\n";

trait Nested
{
    use Counts;
    public function twice()
    {
        return $this->bump()->bump()->count();
    }
}
class UsesNested
{
    use Nested;
}
echo "nested trait: ", (new UsesNested)->twice(), "\n";

class OverridesTrait
{
    use Greets;
    public function greet($who)
    {
        return "class method wins for " . $who;
    }
}
echo (new OverridesTrait)->greet("x"), "\n";

class Parent2
{
    public function hi()
    {
        return "parent hi";
    }
}
trait HiTrait
{
    public function hi()
    {
        return "trait hi over " . parent::hi();
    }
}
class Child2 extends Parent2
{
    use HiTrait;
}
echo (new Child2)->hi(), "\n";
echo "instanceof trait name: ", ($p instanceof Greets ? "yes" : "no"), "\n";
