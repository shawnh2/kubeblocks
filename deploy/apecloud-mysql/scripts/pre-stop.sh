#!/bin/bash
drop_followers() {
echo "leader=$leader" >> /data/mysql/.kb_pre_stop.log
echo "KB_POD_NAME=$KB_POD_NAME" >> /data/mysql/.kb_pre_stop.log
if [ -z "$leader" -o "$KB_POD_NAME" = "$leader" ]; then
  echo "no leader or self is leader, exit" >> /data/mysql/.kb_pre_stop.log
  exit 0
fi
host=$(eval echo \$KB_"$idx"_HOSTNAME)
echo "host=$host" >> /data/mysql/.kb_pre_stop.log
leader_idx=${leader##*-}
leader_host=$(eval echo \$KB_"$leader_idx"_HOSTNAME)
if [ ! -z $leader_host ]; then 
  host_flag="-h$leader_host"
fi
if [ ! -z $MYSQL_ROOT_PASSWORD ]; then 
  password_flag="-p$MYSQL_ROOT_PASSWORD"
fi
echo "mysql $host_flag -uroot $password_flag -e \"call dbms_consensus.downgrade_follower('$host:13306');\" 2>&1 " >> /data/mysql/.kb_pre_stop.log
mysql $host_flag -uroot $password_flag -e "call dbms_consensus.downgrade_follower('$host:13306');" 2>&1
echo "mysql $host_flag -uroot $password_flag -e \"call dbms_consensus.drop_learner('$host:13306');\" 2>&1 " >> /data/mysql/.kb_pre_stop.log
mysql $host_flag -uroot $password_flag -e "call dbms_consensus.drop_learner('$host:13306');" 2>&1
}

set_my_weight_to_zero() {
  if [ ! -z $MYSQL_ROOT_PASSWORD ]; then 
    password_flag="-p$MYSQL_ROOT_PASSWORD"
  fi

  if [ "$KB_POD_NAME" = "$leader" ]; then
    echo "self is leader, before scale in, need to set my election weight to 0." >> /data/mysql/.kb_pre_stop.log
    host=$(eval echo \$KB_"$idx"_HOSTNAME)
    echo "set weight to 0. mysql -uroot $password_flag -e \"call dbms_consensus.configure_follower('$host:13306',0 ,false);\" 2>&1" >> /data/mysql/.kb_pre_stop.log
    mysql -uroot $password_flag -e "call dbms_consensus.configure_follower('$host:13306',0 ,false);" 2>&1
  fi
}

switchover() {
  if [ ! -z $MYSQL_ROOT_PASSWORD ]; then 
    password_flag="-p$MYSQL_ROOT_PASSWORD"
  fi
  #new_leader_host=$KB_0_HOSTNAME
  if [ "$KB_POD_NAME" = "$leader" ]; then
    echo "self is leader, need to switchover" >> /data/mysql/.kb_pre_stop.log
    echo "try to get global cluster info" >> /data/mysql/.kb_pre_stop.log
    global_info=`mysql -uroot $password_flag 2>/dev/null -e "select IP_PORT from information_schema.wesql_cluster_global order by MATCH_INDEX desc;"`
    echo "all nodes: $global_info" >> /data/mysql/.kb_pre_stop.log
    global_info_arr=($global_info)
    echo "all nodes array: ${global_info_arr[0]},${global_info_arr[1]},${global_info_arr[2]},${global_info_arr[3]}"  >> /data/mysql/.kb_pre_stop.log
    echo "array size: ${#global_info_arr[@]}, the first one is not real address,just the field name IP_PORT"  >> /data/mysql/.kb_pre_stop.log

    host=$(eval echo \$KB_"$idx"_HOSTNAME)
    host_ip_port=$host:13306
    try_times=10
    for((i=1;i<${#global_info_arr[@]};i++)) do
      if [ "$host_ip_port" == "${global_info_arr[i]}" ];then
        echo "do not transfer to leader, leader:${global_info_arr[i]}"  >> /data/mysql/.kb_pre_stop.log;
      else
        echo "try to transfer to:${global_info_arr[i]}"  >> /data/mysql/.kb_pre_stop.log;
        echo "mysql -uroot $password_flag -e \"call dbms_consensus.change_leader('${global_info_arr[i]}');\" 2>&1" >> /data/mysql/.kb_pre_stop.log
        mysql -uroot $password_flag -e "call dbms_consensus.change_leader('${global_info_arr[i]}');" 2>&1
        sleep 1
        role_info=`mysql -uroot $password_flag 2>/dev/null -e "select ROLE from information_schema.wesql_cluster_local;"`
        role_info_arr=($role_info)
        real_role=${role_info_arr[1]}
        echo "this node's current role info:$real_role"  >> /data/mysql/.kb_pre_stop.log
        if [ "$real_role" == "Follower" ];then
          echo "transfer successfully" >> /data/mysql/.kb_pre_stop.log
          new_leader_host_and_port=${global_info_arr[i]}
          # get rid of port
          new_leader_host=${new_leader_host_and_port%%:*}
          echo "new_leader_host=$new_leader_host" >> /data/mysql/.kb_pre_stop.log
          leader=`echo "$new_leader_host" | cut -d "." -f 1`
          echo "leader_host: $leader"  >> /data/mysql/.kb_pre_stop.log
          idx=${KB_POD_NAME##*-}
          break
        fi
      fi
      ((try_times--))
      if [ $try_times -le 0 ];then
        echo "try too many times" >> /data/mysql/.kb_pre_stop.log
        break
      fi
    done
  fi
}
leader=`cat /etc/annotations/leader`
idx=${KB_POD_NAME##*-}
current_component_replicas=`cat /etc/annotations/component-replicas`
echo "current replicas: $current_component_replicas" >> /data/mysql/.kb_pre_stop.log
if [ ! $idx -lt $current_component_replicas ] && [ $current_component_replicas -ne 0 ]; then 
    # if idx greater than or equal to current_component_replicas means the cluster's scaling in
    # put .restore on pvc for next scaling out, if pvc not deleted
    touch /data/mysql/data/.restore; sync
    # set wegiht to 0 and switch leader before leader scaling in itself
    set_my_weight_to_zero
    switchover
    # only scaling in need to drop followers
    drop_followers
elif [ $current_component_replicas -eq 0 ]; then
    # stop, do nothing.
    echo "stop, do nothing" >> /data/mysql/.kb_pre_stop.log
else 
    # restart, switchover first.
    echo "Also try to switchover just before restart" >> /data/mysql/.kb_pre_stop.log
    switchover
    echo "no need to drop followers" >> /data/mysql/.kb_pre_stop.log
fi
