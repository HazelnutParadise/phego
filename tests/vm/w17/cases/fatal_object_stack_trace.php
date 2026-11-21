<?php
class Repo
{
    public function find($id, $opts = [])
    {
        return $this->load($id, "row");
    }

    private function load($id, $kind)
    {
        return Loader::fetch($id, 2.5, null, true);
    }
}

class Loader
{
    public static function fetch($id, $f, $n, $b)
    {
        return undefined_helper($id);
    }
}

function run()
{
    $r = new Repo;
    return $r->find(42, ["a" => 1]);
}
echo "running\n";
run();
