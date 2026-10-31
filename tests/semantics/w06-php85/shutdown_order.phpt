--TEST--
Request shutdown destroys solely owned globals in reverse order before remaining objects
--FILE--
<?php
class S { public $name; function __construct($n){$this->name=$n;} function __destruct(){ echo "destruct {$this->name}\n"; } }
$first = new S('first-global');
$shared = new S('shared');
$alias = $shared;
$second = new S('second-global');
$holder = [new S('in-array')];
echo "end of script\n";
--EXPECT--
end of script
destruct second-global
destruct first-global
destruct shared
destruct in-array
