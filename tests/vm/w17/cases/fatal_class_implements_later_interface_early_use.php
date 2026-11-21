<?php
echo "start\n";
echo (new Impl)->m(), "\n";
class Impl implements LaterIface
{
    public function m()
    {
        return "impl";
    }
}
interface LaterIface
{
    public function m();
}
