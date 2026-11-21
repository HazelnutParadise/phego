<?php
class Repo
{
    public function find($id): ?Repo
    {
        return $id > 0 ? $this : "missing";
    }
}
$r = new Repo;
echo ($r->find(1) === $r ? "found" : "no"), "\n";
$r->find(0);
