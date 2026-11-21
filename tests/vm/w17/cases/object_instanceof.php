<?php
interface Walks
{
}
interface Runs extends Walks
{
}
class Animal
{
}
class Dog extends Animal implements Runs
{
}
class Cat extends Animal
{
}

function yn($v)
{
    return $v ? "yes" : "no";
}

$d = new Dog;
$c = new Cat;
echo "dog instanceof Dog ", yn($d instanceof Dog), "\n";
echo "dog instanceof Animal ", yn($d instanceof Animal), "\n";
echo "dog instanceof Runs ", yn($d instanceof Runs), "\n";
echo "dog instanceof Walks ", yn($d instanceof Walks), "\n";
echo "cat instanceof Walks ", yn($c instanceof Walks), "\n";
echo "cat instanceof Dog ", yn($c instanceof Dog), "\n";
echo "negated ", yn(!$c instanceof Dog), "\n";

$cls = "Animal";
echo "string variable ", yn($d instanceof $cls), "\n";
$cls = "animal";
echo "string variable lower ", yn($d instanceof $cls), "\n";
$other = new Cat;
echo "object on right ", yn($c instanceof $other), " ", yn($d instanceof $other), "\n";

echo "undefined class on right ", yn($d instanceof NoSuchClass), "\n";
$missing = "AlsoMissing";
echo "undefined class string ", yn($d instanceof $missing), "\n";

$i = 5;
$s = "Dog";
$n = null;
$a = [1];
echo "int left ", yn($i instanceof Dog), "\n";
echo "string left ", yn($s instanceof Dog), "\n";
echo "null left ", yn($n instanceof Dog), "\n";
echo "array left ", yn($a instanceof Dog), "\n";

class Self_
{
    public function check($o)
    {
        return yn($o instanceof self) . yn($o instanceof static);
    }
}
class SelfChild extends Self_
{
}
echo "self/static: ", (new SelfChild)->check(new Self_), " ", (new SelfChild)->check(new SelfChild), "\n";
