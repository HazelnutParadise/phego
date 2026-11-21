<?php
class Vault
{
    public function open()
    {
        return "opened with " . $this->code();
    }

    private function code()
    {
        return "1234";
    }
}
$v = new Vault;
echo $v->open(), "\n";
echo $v->code(), "\n";
