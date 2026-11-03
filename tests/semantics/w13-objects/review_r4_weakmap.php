<?php
// REQ-018/021; T-035/036/041/042.
class W13R4Stringable { public function __toString(): string { $GLOBALS['r4_calls']++; return 'value'; } }
class W13R4Ordinary {}
class W13R4WeakMapLazy { public int $p; }


$w13R4Names = ['empty','sameentries','clone','differentkey','differentvalue','differentcount','reversedinsertorder','arrayvalues','selfcycle','mutualcycle','callback-values','sharedidentity','ordinary-class'];
foreach ($w13R4Names as $case) {
    $tests['r4-weakmap-'.$case] = function() use ($case) {
        $GLOBALS['r4_calls'] = 0;
        $a = new WeakMap; $b = new WeakMap;
        $k1 = new stdClass; $k2 = new stdClass;
        switch ($case) {
            case 'sameentries': $a[$k1]=1; $b[$k1]=1; break;
            case 'clone': $a[$k1]=1; $b=clone $a; break;
            case 'differentkey': $a[$k1]=1; $b[$k2]=1; break;
            case 'differentvalue': $a[$k1]=1; $b[$k1]=2; break;
            case 'differentcount': $a[$k1]=1; $b[$k1]=1; $b[$k2]=2; break;
            case 'reversedinsertorder': $a[$k1]=1; $a[$k2]=2; $b[$k2]=2; $b[$k1]=1; break;
            case 'arrayvalues': $a[$k1]=[1]; $b[$k1]=[2]; break;
            case 'selfcycle': $a[$k1]=$a; $b[$k1]=$b; break;
            case 'mutualcycle': $a[$k1]=$b; $b[$k2]=$a; break;
            case 'callback-values': $a[$k1]=new W13R4Stringable; $b[$k1]=new W13R4Stringable; $r=new ReflectionClass(W13R4WeakMapLazy::class); $a[$k2]=$r->newLazyGhost(function() { $GLOBALS['r4_calls']++; throw new Exception('lazy initializer called'); }); $b[$k2]=$r->newLazyGhost(function() { $GLOBALS['r4_calls']++; throw new Exception('lazy initializer called'); }); break;
            case 'sharedidentity': $b=$a; break;
            case 'ordinary-class': $b=new W13R4Ordinary; break;
        }
        return [attempt(fn() => $a <=> $b), attempt(fn() => $b <=> $a), attempt(fn() => $a == $b), attempt(fn() => $a === $b), $GLOBALS['r4_calls']];
    };
}
