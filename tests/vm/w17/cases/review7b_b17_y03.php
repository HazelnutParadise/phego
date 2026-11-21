<?php
trait T { abstract function m(self $x): self; }
class C { use T; function m(self $x): self { return $this; } }
echo "ok\n";
