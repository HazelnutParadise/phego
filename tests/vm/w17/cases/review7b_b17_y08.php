<?php
class P {}
trait T { abstract function m(parent $x); }
class C extends P { use T; function m(P $x) {} }
echo "ok\n";
