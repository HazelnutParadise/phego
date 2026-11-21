<?php
trait T { function m() {} }
class A { use T { Nope::zz as yy; } }
echo "x\n";
