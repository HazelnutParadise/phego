<?php
echo "start\n";
class Quiet
{
    private function __call($n, $a)
    {
        return "private __call still used for " . $n;
    }

    protected function __get($n)
    {
        return "protected __get still used for " . $n;
    }

    private static function __callStatic($n, $a)
    {
        return "private __callStatic still used for " . $n;
    }
}
$q = new Quiet;
echo $q->anything(), "\n";
echo $q->prop, "\n";
echo Quiet::stat(), "\n";
