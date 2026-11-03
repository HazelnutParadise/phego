<?php
// REQ-018/020; T-035/036/039/040. Initialized proxies retain wrapper hook dispatch.
class W13R10HookParent {
    public int $spare = 2;
    public int $x = 1 {
        &get { if (false) return $this->x; $GLOBALS['w13_r10_hook_events'][] = 'parent:get'; return $GLOBALS['w13_r10_parent_cell']; }
    }
    public int $y {
        set { $GLOBALS['w13_r10_hook_events'][] = ['parent:set', $value]; }
    }
}
class W13R10HookChild extends W13R10HookParent {
    public int $x = 1 {
        &get { if (false) return $this->x; $GLOBALS['w13_r10_hook_events'][] = 'wrapper:get'; return $GLOBALS['w13_r10_child_cell']; }
    }
    public int $y {
        set { $GLOBALS['w13_r10_hook_events'][] = ['wrapper:set', $value]; }
    }
}
$w13R10ProxyHookNames = [];
foreach ([false, true] as $warm) foreach (['read', 'write', 'reference', 'isset'] as $operation) {
    $name = 'r10-proxy-hooks-'.($warm ? 'warm' : 'cold').'-'.$operation;
    $w13R10ProxyHookNames[] = $name;
    $tests[$name] = function() use($warm, $operation) {
        $GLOBALS['w13_r10_hook_events'] = [];
        $GLOBALS['w13_r10_parent_cell'] = 11; $GLOBALS['w13_r10_child_cell'] = 7;
        $backing = new W13R10HookParent; $reflection = new ReflectionClass(W13R10HookChild::class);
        $proxy = $reflection->newLazyProxy(function() use($backing) { $GLOBALS['w13_r10_hook_events'][] = 'factory'; return $backing; });
        if ($warm) $GLOBALS['w13_r10_hook_events'][] = ['warm', $proxy->spare];
        $result = w13R10Attempt(function() use($proxy, $operation) {
            switch ($operation) {
                case 'read': return $proxy->x;
                case 'write': $proxy->y = 3; return null;
                case 'reference': $alias =& $proxy->x; $alias = 3; return $alias;
                case 'isset': return isset($proxy->x);
            }
        });
        return [$result, $GLOBALS['w13_r10_child_cell'], $GLOBALS['w13_r10_parent_cell'], $GLOBALS['w13_r10_hook_events'], $reflection->isUninitializedLazyObject($proxy)];
    };
}
