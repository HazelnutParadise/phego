<?php
class A
{
    public function make(): static
    {
        return new A;
    }
}
class B extends A
{
}
echo (new A)->make()::class, "\n";
(new B)->make();
