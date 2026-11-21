<?php
class Bad
{
    public function __destruct()
    {
        echo "destructor runs\n";
        $this->missing();
    }
}
$b = new Bad;
echo "unsetting\n";
unset($b);
echo "not reached\n";
