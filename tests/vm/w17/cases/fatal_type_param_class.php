<?php
class Cat
{
}
class Dog
{
}
class Vet
{
    public function treat(Dog $d)
    {
        return "treated";
    }
}
$v = new Vet;
echo $v->treat(new Dog), "\n";
echo $v->treat(new Cat), "\n";
