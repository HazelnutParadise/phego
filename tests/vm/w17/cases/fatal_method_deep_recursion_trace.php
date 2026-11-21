<?php
class Walker
{
    public function walk($n)
    {
        if ($n === 0) {
            return $this->missing($n);
        }
        return $this->walk($n - 1);
    }
}
echo "walking\n";
(new Walker)->walk(3);
