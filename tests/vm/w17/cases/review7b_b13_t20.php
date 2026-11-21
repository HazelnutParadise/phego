<?php
trait T { abstract function m(self $a); }
class C { use T; function m(C $a) {} }
echo "ok\n";
