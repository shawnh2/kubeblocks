/*
Copyright ApeCloud Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dataprotection

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sethvargo/go-password/password"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/util/yaml"

	dataprotectionv1alpha1 "github.com/apecloud/kubeblocks/apis/dataprotection/v1alpha1"
	intctrlutil "github.com/apecloud/kubeblocks/internal/controllerutil"
	testdbaas "github.com/apecloud/kubeblocks/internal/testutil/dbaas"
)

var _ = Describe("BackupPolicyTemplate Controller", func() {

	const waitDuration = time.Second * 3

	var ctx = context.Background()

	cleanEnv := func() {
		// must wait until resources deleted and no longer exist before the testcases start,
		// otherwise if later it needs to create some new resource objects with the same name,
		// in race conditions, it will find the existence of old objects, resulting failure to
		// create the new objects.
		By("clean resources")

		// delete rest mocked objects
		ml := client.HasLabels{testCtx.TestObjLabelKey}

		// non-namespaced
		testdbaas.ClearResources(&testCtx, intctrlutil.BackupToolSignature, ml)
		testdbaas.ClearResources(&testCtx, intctrlutil.BackupPolicyTemplateSignature, ml)
	}

	BeforeEach(cleanEnv)

	AfterEach(cleanEnv)

	AfterEach(func() {
		// Add any teardown steps that needs to be executed after each test
	})

	genarateNS := func(prefix string) types.NamespacedName {
		randomStr, _ := password.Generate(6, 0, 0, true, false)
		key := types.NamespacedName{
			Name:      prefix + randomStr,
			Namespace: "default",
		}
		return key
	}

	assureBackupPolicyTemplateObj := func(backupTool string) *dataprotectionv1alpha1.BackupPolicyTemplate {
		By("By assure an backupPolicyTemplate obj")
		backupPolicyYaml := `
apiVersion: dataprotection.kubeblocks.io/v1alpha1
kind: BackupPolicyTemplate
metadata:
  name: backup-policy-template-demo
  namespace: default
spec:
  schedule: "0 3 * * *"
  ttl: 168h0m0s
  backupToolName: xtrabackup-mysql
  onFailAttempted: 3
`
		backupPolicyTemp := &dataprotectionv1alpha1.BackupPolicyTemplate{}
		Expect(yaml.Unmarshal([]byte(backupPolicyYaml), backupPolicyTemp)).Should(Succeed())
		ns := genarateNS("backup-policy-template-")
		backupPolicyTemp.Name = ns.Name
		backupPolicyTemp.Namespace = ns.Namespace
		backupPolicyTemp.Spec.BackupToolName = backupTool
		Expect(testCtx.CreateObj(ctx, backupPolicyTemp)).Should(Succeed())
		return backupPolicyTemp
	}

	assureBackupToolObj := func() *dataprotectionv1alpha1.BackupTool {
		By("By assure an backupTool obj")
		backupToolYaml := `
apiVersion: dataprotection.kubeblocks.io/v1alpha1
kind: BackupTool
metadata:
  name: xtrabackup-mysql
  namespace: default
spec:
  image: percona/percona-xtrabackup
  databaseEngine: mysql
  deployKind: job
  resources:
    limits:
      cpu: "1"
      memory: 2Gi

  env:
    - name: DATA_DIR
      value: /var/lib/mysql

    - name: BACKUP_DIR_PREFIX
      valueFrom:
        fieldRef:
          fieldPath: metadata.namespace

    - name: BACKUP_DIR
      value: /data/$(BACKUP_DIR_PREFIX)

  restoreCommands:
    - |
      echo "BACKUP_DIR=${BACKUP_DIR} BACKUP_NAME=${BACKUP_NAME} DATA_DIR=${DATA_DIR}" && \
      mkdir -p /tmp/data/ && cd /tmp/data \
      && xbstream -x < /${BACKUP_DIR}/${BACKUP_NAME}.xbstream \
      && xtrabackup --decompress  --target-dir=/tmp/data/ \
      && find . -name "*.qp"|xargs rm -f \
      && rm -rf ${DATA_DIR}/* \
      && rsync -avrP /tmp/data/ ${DATA_DIR}/ \
      && rm -rf /tmp/data/ \
      && chmod -R 0777 ${DATA_DIR}

  # Optional
  incrementalRestoreCommands: []
  backupCommands:
    - echo "DB_HOST=${DB_HOST} DB_USER=${DB_USER} DB_PASSWORD=${DB_PASSWORD} DATA_DIR=${DATA_DIR} BACKUP_DIR=${BACKUP_DIR} BACKUP_NAME=${BACKUP_NAME}";
      mkdir -p /${BACKUP_DIR};
      xtrabackup --compress --backup  --safe-slave-backup --slave-info --stream=xbstream --host=${DB_HOST} --user=${DB_USER} --password=${DB_PASSWORD} --datadir=${DATA_DIR} > /${BACKUP_DIR}/${BACKUP_NAME}.xbstream
  # Optional
  incrementalBackupCommands: []
`
		backupTool := &dataprotectionv1alpha1.BackupTool{}
		Expect(yaml.Unmarshal([]byte(backupToolYaml), backupTool)).Should(Succeed())
		ns := genarateNS("backup-tool-")
		backupTool.Name = ns.Name
		backupTool.Namespace = ns.Namespace
		Expect(testCtx.CreateObj(ctx, backupTool)).Should(Succeed())
		return backupTool
	}

	Context("When creating backupPolicyTemplate", func() {
		It("Should success with no error", func() {

			By("By creating a backupTool")
			backupTool := assureBackupToolObj()

			By("By creating a backupPolicyTemplate from backupTool: " + backupTool.Name)
			toCreate := assureBackupPolicyTemplateObj(backupTool.Name)
			key := types.NamespacedName{
				Name:      toCreate.Name,
				Namespace: toCreate.Namespace,
			}
			time.Sleep(waitDuration)
			result := &dataprotectionv1alpha1.BackupPolicyTemplate{}
			Expect(k8sClient.Get(ctx, key, result)).Should(Succeed())
		})
	})

})
