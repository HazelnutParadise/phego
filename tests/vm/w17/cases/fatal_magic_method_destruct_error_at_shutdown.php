<?php
class Bad
{
    public function __destruct()
    {
        echo "shutdown destructor\n";
        echo 1 % 0;
    }
}
class Good
{
    public function __destruct()
    {
        echo "good destructor\n";
    }
}
$g = new Good;
$b = new Bad;
echo "end of script\n";
