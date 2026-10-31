--TEST--
Destructors that release children run nested
--FILE--
<?php
class E { public $child; public $name; function __construct($n,$c=null){$this->name=$n;$this->child=$c;} function __destruct(){ echo "enter {$this->name}\n"; $this->child = null; echo "leave {$this->name}\n"; } }
$top = new E('top', new E('mid', new E('leaf')));
unset($top);
echo "done\n";
--EXPECT--
enter top
enter mid
enter leaf
leave leaf
leave mid
leave top
done
