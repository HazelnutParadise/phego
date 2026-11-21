<?php
class OnlyInstance
{
    public function __call($n, $a)
    {
        return "instance " . $n;
    }
}
echo (new OnlyInstance)->thing(), "\n";
echo OnlyInstance::thing(), "\n";
