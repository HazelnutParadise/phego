<?php
function log_it($m): void
{
    echo "log: ", $m, "\n";
    if ($m === "early") {
        return;
    }
    echo "after\n";
}
$r = log_it("first");
echo "void returns null: ", ($r === null ? "yes" : "no"), "\n";
log_it("early");

class Svc
{
    public function run(): void
    {
        echo "run\n";
    }
    public function stop(): never
    {
        echo "stopping\n";
        exit(3);
    }
}
(new Svc)->run();
echo "before never\n";
(new Svc)->stop();
echo "not reached\n";
