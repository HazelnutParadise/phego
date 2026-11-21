<?php
trait T { abstract function m(): self; }
class C { use T; function m(): C { return $this; } }
echo "ok\n";
