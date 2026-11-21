<?php
echo "start\n";
trait NeedsLabel
{
    abstract public function label();
    public function show()
    {
        return $this->label();
    }
}
class NoLabel
{
    use NeedsLabel;
}
